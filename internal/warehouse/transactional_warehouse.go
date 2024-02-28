package warehouse

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kiwicom/terraform-provider-montecarlo/client"
	"github.com/kiwicom/terraform-provider-montecarlo/internal/common"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &TransactionalWarehouseResource{}
var _ resource.ResourceWithImportState = &TransactionalWarehouseResource{}

// To simplify provider implementations, a named function can be created with the resource implementation.
func NewTransactionalWarehouseResource() resource.Resource {
	return &TransactionalWarehouseResource{}
}

// TransactionalWarehouseResource defines the resource implementation.
type TransactionalWarehouseResource struct {
	client client.MonteCarloClient
}

// TransactionalWarehouseResourceModel describes the resource data model according to its Schema.
type TransactionalWarehouseResourceModel struct {
	Uuid               types.String             `tfsdk:"uuid"`
	Name               types.String             `tfsdk:"name"`
	DbType             types.String             `tfsdk:"db_type"`
	CollectorUuid      types.String             `tfsdk:"collector_uuid"`
	Credentials        TransactionalCredentials `tfsdk:"credentials"`
	DeletionProtection types.Bool               `tfsdk:"deletion_protection"`
}

type TransactionalCredentials struct {
	ConnectionUuid types.String `tfsdk:"connection_uuid"`
	Host           types.String `tfsdk:"host"`
	Port           types.Int64  `tfsdk:"port"`
	Database       types.String `tfsdk:"database"`
	Username       types.String `tfsdk:"username"`
	Password       types.String `tfsdk:"password"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (m TransactionalWarehouseResourceModel) GetUuid() types.String          { return m.Uuid }
func (m TransactionalWarehouseResourceModel) GetCollectorUuid() types.String { return m.CollectorUuid }
func (m TransactionalWarehouseResourceModel) GetName() types.String          { return m.Name }
func (m TransactionalWarehouseResourceModel) GetConnectionUuid() types.String {
	return m.Credentials.ConnectionUuid
}

type TransactionalWarehouseResourceModelVO struct {
	Uuid               types.String    `tfsdk:"uuid"`
	ConnectionUuid     types.String    `tfsdk:"connection_uuid"`
	Name               types.String    `tfsdk:"name"`
	DbType             types.String    `tfsdk:"db_type"`
	CollectorUuid      types.String    `tfsdk:"collector_uuid"`
	Configuration      ConfigurationV0 `tfsdk:"configuration"`
	DeletionProtection types.Bool      `tfsdk:"deletion_protection"`
}

type ConfigurationV0 struct {
	Host     types.String `tfsdk:"host"`
	Port     types.Int64  `tfsdk:"port"`
	Database types.String `tfsdk:"database"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

func (r *TransactionalWarehouseResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_transactional_warehouse"
}

func (r *TransactionalWarehouseResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		Attributes: map[string]schema.Attribute{
			"uuid": schema.StringAttribute{
				Computed: true,
				Optional: false,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"db_type": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf("POSTGRES", "MYSQL", "SQL-SERVER"),
				},
			},
			"collector_uuid": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"credentials": schema.SingleNestedAttribute{
				Required: true,
				Attributes: map[string]schema.Attribute{
					"connection_uuid": schema.StringAttribute{
						Computed: true,
						Optional: false,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"host": schema.StringAttribute{
						Required: true,
					},
					"port": schema.Int64Attribute{
						Required: true,
					},
					"database": schema.StringAttribute{
						Required: true,
					},
					"username": schema.StringAttribute{
						Required:  true,
						Sensitive: true,
					},
					"password": schema.StringAttribute{
						Required:  true,
						Sensitive: true,
					},
					"updated_at": schema.StringAttribute{
						Computed: true,
						Optional: false,
					},
				},
			},
			"deletion_protection": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
		},
	}
}

func (r *TransactionalWarehouseResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := common.Configure(req)
	resp.Diagnostics.Append(diags...)
	r.client = client
}

func (r *TransactionalWarehouseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TransactionalWarehouseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, diags := addConnection(ctx, r.client, r, data, client.TrxConnectionType, TrxKeyExtractor)
	resp.Diagnostics.Append(diags...)
	if result == nil {
		return
	}

	data.Uuid = types.StringValue(result.AddConnection.Connection.Warehouse.Uuid)
	data.Credentials.UpdatedAt = types.StringValue(result.AddConnection.Connection.CreatedOn)
	data.Credentials.ConnectionUuid = types.StringValue(result.AddConnection.Connection.Uuid)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TransactionalWarehouseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TransactionalWarehouseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	getResult := client.GetWarehouse{}
	variables := map[string]interface{}{"uuid": client.UUID(data.Uuid.ValueString())}

	if bytes, err := r.client.ExecRaw(ctx, client.GetWarehouseQuery, variables); err != nil && len(bytes) == 0 {
		toPrint := fmt.Sprintf("MC client 'GetWarehouse' query result - %s", err.Error())
		resp.Diagnostics.AddError(toPrint, "")
		return
	} else if jsonErr := json.Unmarshal(bytes, &getResult); jsonErr != nil {
		toPrint := fmt.Sprintf("MC client 'GetWarehouse' query failed to unmarshal data - %s", jsonErr.Error())
		resp.Diagnostics.AddError(toPrint, "")
		return
	} else if getResult.GetWarehouse == nil {
		toPrint := fmt.Sprintf("MC client 'GetWarehouse' query failed to find warehouse [uuid: %s]. "+
			"This resource will be removed from the Terraform state without deletion.", data.Uuid.ValueString())
		if err != nil {
			toPrint = fmt.Sprintf("%s - %s", toPrint, err.Error())
		} // response missing warehouse data may or may not contain error
		resp.Diagnostics.AddWarning(toPrint, "")
		resp.State.RemoveResource(ctx)
		return
	}

	readCollectorUuid := getResult.GetWarehouse.DataCollector.Uuid
	confCollectorUuid := data.CollectorUuid.ValueString()
	if readCollectorUuid != confCollectorUuid {
		resp.Diagnostics.AddWarning(fmt.Sprintf("Obtained Transactional warehouse with [uuid: %s] but its Data "+
			"Collector UUID does not match with configured value [obtained: %s, configured: %s]. Transactional "+
			"warehouse might have been moved to other Data Collector externally. This resource will be removed "+
			"from the Terraform state without deletion.",
			data.Uuid.ValueString(), readCollectorUuid, confCollectorUuid), "")
		resp.State.RemoveResource(ctx)
		return
	}

	readCredentials := TransactionalCredentials{
		ConnectionUuid: types.StringNull(),
		Host:           types.StringNull(),
		Port:           types.Int64Null(),
		Database:       types.StringNull(),
		Username:       types.StringNull(),
		Password:       types.StringNull(),
		UpdatedAt:      types.StringNull(),
	}

	for _, connection := range getResult.GetWarehouse.Connections {
		if connection.Uuid == data.Credentials.ConnectionUuid.ValueString() {
			if connection.Type != client.TrxConnectionTypeResponse {
				resp.Diagnostics.AddError(
					fmt.Sprintf("Obtained Warehouse [uuid: %s, connection_uuid: %s] but got unexpected connection "+
						"type '%s'", data.Uuid.ValueString(), connection.Uuid, connection.Type),
					"Users can manually fix remote state or delete this resource from the Terraform configuration.")
				return
			}

			readCredentials = data.Credentials
			readCredentials.UpdatedAt = types.StringValue(connection.UpdatedOn)
			if connection.UpdatedOn == "" {
				readCredentials.UpdatedAt = types.StringValue(connection.CreatedOn)
			}
		}
	}

	if !readCredentials.ConnectionUuid.IsNull() && !readCredentials.UpdatedAt.Equal(data.Credentials.UpdatedAt) {
		readCredentials.Host = types.StringValue("(unknown remote value)")
		readCredentials.Port = types.Int64Value(-1)
		readCredentials.Database = types.StringValue("(unknown remote value)")
		readCredentials.Username = types.StringValue("(unknown remote value)")
		readCredentials.Password = types.StringValue("(unknown remote value)")
	}

	data.Credentials = readCredentials
	data.Name = types.StringValue(getResult.GetWarehouse.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TransactionalWarehouseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TransactionalWarehouseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	setNameResult := client.SetWarehouseName{}
	variables := map[string]interface{}{
		"dwId": client.UUID(data.Uuid.ValueString()),
		"name": data.Name.ValueString(),
	}

	if err := r.client.Mutate(ctx, &setNameResult, variables); err != nil {
		to_print := fmt.Sprintf("MC client 'SetWarehouseName' mutation result - %s", err.Error())
		resp.Diagnostics.AddError(to_print, "")
		return
	}

	if data.Credentials.ConnectionUuid.IsUnknown() || data.Credentials.ConnectionUuid.IsNull() {
		if result, diags := addConnection(ctx, r.client, r, data, client.TrxConnectionType, TrxKeyExtractor); result != nil {
			resp.Diagnostics.Append(diags...)
			data.Credentials.UpdatedAt = types.StringValue(result.AddConnection.Connection.CreatedOn)
			data.Credentials.ConnectionUuid = types.StringValue(result.AddConnection.Connection.Uuid)
		} else {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	if updateResult, diags := updateConnection(ctx, r.client, r, data, TrxKeyExtractor); updateResult == nil {
		resp.Diagnostics.Append(diags...)
	} else {
		data.Credentials.UpdatedAt = types.StringValue(updateResult.UpdateCredentialsV2.UpdatedAt)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	}
}

func (r *TransactionalWarehouseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TransactionalWarehouseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.DeletionProtection.ValueBool() {
		resp.Diagnostics.AddError(
			"Failed to delete warehouse because deletion_protection is set to true. "+
				"Set it to false to proceed with warehouse deletion",
			"Deletion protection flag will prevent this resource deletion even if it was already deleted "+
				"from the real system. For reasons why this is preferred behaviour check out documentation.",
		)
		return
	}

	removeResult := client.RemoveConnection{}
	variables := map[string]interface{}{"connectionId": client.UUID(data.Credentials.ConnectionUuid.ValueString())}
	if err := r.client.Mutate(ctx, &removeResult, variables); err != nil {
		toPrint := fmt.Sprintf("MC client 'RemoveConnection' mutation result - %s", err.Error())
		resp.Diagnostics.AddError(toPrint, "")
	} else if !removeResult.RemoveConnection.Success {
		toPrint := "MC client 'RemoveConnection' mutation - success = false, " +
			"connection probably already doesn't exists. This resource will continue with its deletion"
		resp.Diagnostics.AddWarning(toPrint, "")
	}
}

func (r *TransactionalWarehouseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idsImported := strings.Split(req.ID, ",")
	if len(idsImported) == 3 && idsImported[0] != "" && idsImported[1] != "" && idsImported[2] != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), idsImported[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("credentials").AtName("connection_uuid"), idsImported[1])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("collector_uuid"), idsImported[2])...)
	} else {
		resp.Diagnostics.AddError("Unexpected Import Identifier", fmt.Sprintf(
			"Expected import identifier with format: <warehouse_uuid>,<connection_uuid>,<data_collector_uuid>. Got: %q", req.ID),
		)
	}
}

func (r *TransactionalWarehouseResource) testCredentials(ctx context.Context, data TransactionalWarehouseResourceModel) (*client.TestDatabaseCredentials, diag.Diagnostics) {
	var diagsResult diag.Diagnostics
	testResult := client.TestDatabaseCredentials{}
	variables := map[string]interface{}{
		"connectionType": client.TrxConnectionType,
		"dbType":         strings.ToLower(data.DbType.ValueString()),
		"host":           data.Credentials.Host.ValueString(),
		"port":           data.Credentials.Port.ValueInt64(),
		"dbName":         data.Credentials.Database.ValueString(),
		"user":           data.Credentials.Username.ValueString(),
		"password":       data.Credentials.Password.ValueString(),
	}

	if err := r.client.Mutate(ctx, &testResult, variables); err != nil {
		toPrint := fmt.Sprintf("MC client 'TestDatabaseCredentials' mutation result - %s", err.Error())
		diagsResult.AddError(toPrint, "")
		return nil, diagsResult
	} else if !testResult.TestDatabaseCredentials.Success {
		diags := databaseTestDiagnosticsToDiags(testResult.TestDatabaseCredentials.Warnings)
		diags = append(diags, databaseTestDiagnosticsToDiags(testResult.TestDatabaseCredentials.Validations)...)
		diagsResult.Append(diags...)
		return nil, diagsResult
	} else {
		return &testResult, diagsResult
	}
}

func databaseTestDiagnosticsToDiags(in []client.DatabaseTestDiagnostic) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, value := range in {
		diags.AddWarning(value.Message, value.Type)
	}
	return diags
}

func (r *TransactionalWarehouseResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"uuid": schema.StringAttribute{
						Computed: true,
						Optional: false,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"connection_uuid": schema.StringAttribute{
						Computed: true,
						Optional: false,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"name": schema.StringAttribute{
						Required: true,
					},
					"db_type": schema.StringAttribute{
						Required: true,
						Validators: []validator.String{
							stringvalidator.OneOf("POSTGRES", "MYSQL", "SQL-SERVER"),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplaceIfConfigured(),
						},
					},
					"collector_uuid": schema.StringAttribute{
						Required: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplaceIfConfigured(),
						},
					},
					"configuration": schema.SingleNestedAttribute{
						Required: true,
						Attributes: map[string]schema.Attribute{
							"host": schema.StringAttribute{
								Required: true,
							},
							"port": schema.Int64Attribute{
								Required: true,
							},
							"database": schema.StringAttribute{
								Required: true,
								PlanModifiers: []planmodifier.String{
									stringplanmodifier.RequiresReplaceIfConfigured(),
								},
							},
							"username": schema.StringAttribute{
								Required:  true,
								Sensitive: true,
							},
							"password": schema.StringAttribute{
								Required:  true,
								Sensitive: true,
							},
						},
					},
					"deletion_protection": schema.BoolAttribute{
						Optional: true,
						Computed: true,
						Default:  booldefault.StaticBool(true),
					},
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var priorStateData TransactionalWarehouseResourceModelVO
				resp.Diagnostics.Append(req.State.Get(ctx, &priorStateData)...)
				if !resp.Diagnostics.HasError() {
					upgradedStateData := TransactionalWarehouseResourceModel{
						Uuid:          priorStateData.Uuid,
						CollectorUuid: priorStateData.CollectorUuid,
						Name:          priorStateData.Name,
						DbType:        priorStateData.DbType,
						Credentials: TransactionalCredentials{
							ConnectionUuid: priorStateData.ConnectionUuid,
							Host:           priorStateData.Configuration.Host,
							Port:           priorStateData.Configuration.Port,
							Database:       priorStateData.Configuration.Database,
							Username:       priorStateData.Configuration.Username,
							Password:       priorStateData.Configuration.Password,
							UpdatedAt:      types.StringNull(),
						},
					}
					resp.Diagnostics.Append(resp.State.Set(ctx, upgradedStateData)...)
				}
			},
		},
	}
}

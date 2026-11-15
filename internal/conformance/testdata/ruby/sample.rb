require "json"

# CONF_LINE_COMMENT

# CONF_DOC_COMMENT
class ConfWidget
  include Comparable
  attr_reader :conf_name

  def conf_public
    a = "CONF_BODY_A"
    a
  end

  def conf_splat(*conf_args)
    1
  end

  private

  # CONF_PRIVATE_DOC
  def conf_private
    JSON.generate([])
  end

  public

  def conf_other
    b = "CONF_BODY_B"
    b
  end
end
